package federation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

// testClientProvider implements ClientProvider for testing.
type testClientProvider struct {
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	restConfig    *rest.Config
	err           error
}

func (p *testClientProvider) GetClientsForUser(_ context.Context, _ *UserInfo) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	if p.err != nil {
		return nil, nil, nil, p.err
	}
	return p.clientset, p.dynamicClient, p.restConfig, nil
}

func TestValidateAccessCheck(t *testing.T) {
	tests := []struct {
		name    string
		check   *AccessCheck
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil check",
			check:   nil,
			wantErr: true,
			errMsg:  "check is nil",
		},
		{
			name:    "empty verb",
			check:   &AccessCheck{Resource: "pods"},
			wantErr: true,
			errMsg:  "verb is required",
		},
		{
			name:    "empty resource",
			check:   &AccessCheck{Verb: "get"},
			wantErr: true,
			errMsg:  "resource is required",
		},
		{
			name:    "invalid verb",
			check:   &AccessCheck{Verb: "invalid", Resource: "pods"},
			wantErr: true,
			errMsg:  "unknown verb",
		},
		{
			name:    "valid get pods",
			check:   &AccessCheck{Verb: "get", Resource: "pods"},
			wantErr: false,
		},
		{
			name:    "valid list deployments",
			check:   &AccessCheck{Verb: "list", Resource: "deployments", APIGroup: "apps"},
			wantErr: false,
		},
		{
			name:    "valid delete with namespace",
			check:   &AccessCheck{Verb: "delete", Resource: "pods", Namespace: "default"},
			wantErr: false,
		},
		{
			name:    "valid create with all fields",
			check:   &AccessCheck{Verb: "create", Resource: "pods", Namespace: "default", Name: "my-pod"},
			wantErr: false,
		},
		{
			name:    "valid wildcard verb",
			check:   &AccessCheck{Verb: "*", Resource: "pods"},
			wantErr: false,
		},
		{
			name:    "valid watch",
			check:   &AccessCheck{Verb: "watch", Resource: "pods"},
			wantErr: false,
		},
		{
			name:    "valid patch",
			check:   &AccessCheck{Verb: "patch", Resource: "deployments", APIGroup: "apps"},
			wantErr: false,
		},
		{
			name:    "valid deletecollection",
			check:   &AccessCheck{Verb: "deletecollection", Resource: "pods"},
			wantErr: false,
		},
		{
			name:    "valid impersonate",
			check:   &AccessCheck{Verb: "impersonate", Resource: "users"},
			wantErr: false,
		},
		{
			name:    "valid subresource",
			check:   &AccessCheck{Verb: "create", Resource: "pods", Subresource: "exec"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccessCheck(tt.check)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAccessCheck() expected error but got nil")
					return
				}
				if !errors.Is(err, ErrInvalidAccessCheck) {
					t.Errorf("ValidateAccessCheck() error = %v, want ErrInvalidAccessCheck", err)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateAccessCheck() error = %v, want to contain %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAccessCheck() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestManager_CheckAccess(t *testing.T) {
	validUser := &UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}

	tests := []struct {
		name          string
		clusterName   string
		user          *UserInfo
		check         *AccessCheck
		setupClient   func(*fake.Clientset)
		wantAllowed   bool
		wantDenied    bool
		wantReason    string
		wantErr       bool
		wantErrIs     error
		providerErr   error
		closedManager bool
	}{
		{
			name:        "allowed - get pods",
			clusterName: "",
			user:        validUser,
			check:       &AccessCheck{Verb: "get", Resource: "pods", Namespace: "default"},
			setupClient: func(cs *fake.Clientset) {
				cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: true,
							Reason:  "RBAC: allowed by ClusterRoleBinding",
						},
					}, nil
				})
			},
			wantAllowed: true,
			wantReason:  "RBAC: allowed by ClusterRoleBinding",
			wantErr:     false,
		},
		{
			name:        "denied - delete pods",
			clusterName: "",
			user:        validUser,
			check:       &AccessCheck{Verb: "delete", Resource: "pods", Namespace: "production"},
			setupClient: func(cs *fake.Clientset) {
				cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: false,
							Denied:  true,
							Reason:  "RBAC: delete denied",
						},
					}, nil
				})
			},
			wantAllowed: false,
			wantDenied:  true,
			wantReason:  "RBAC: delete denied",
			wantErr:     false,
		},
		{
			name:        "nil user",
			clusterName: "",
			user:        nil,
			check:       &AccessCheck{Verb: "get", Resource: "pods"},
			wantErr:     true,
			wantErrIs:   ErrUserInfoRequired,
		},
		{
			name:        "invalid check - nil",
			clusterName: "",
			user:        validUser,
			check:       nil,
			wantErr:     true,
			wantErrIs:   ErrInvalidAccessCheck,
		},
		{
			name:        "invalid check - empty verb",
			clusterName: "",
			user:        validUser,
			check:       &AccessCheck{Verb: "", Resource: "pods"},
			wantErr:     true,
			wantErrIs:   ErrInvalidAccessCheck,
		},
		{
			name:        "invalid cluster name",
			clusterName: "../../../etc/passwd",
			user:        validUser,
			check:       &AccessCheck{Verb: "get", Resource: "pods"},
			wantErr:     true,
			wantErrIs:   ErrInvalidClusterName,
		},
		{
			name:        "client provider error",
			clusterName: "",
			user:        validUser,
			check:       &AccessCheck{Verb: "get", Resource: "pods"},
			providerErr: errors.New("connection refused"),
			wantErr:     true,
			wantErrIs:   ErrAccessCheckFailed,
		},
		{
			name:        "API server error",
			clusterName: "",
			user:        validUser,
			check:       &AccessCheck{Verb: "get", Resource: "pods"},
			setupClient: func(cs *fake.Clientset) {
				cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("internal server error")
				})
			},
			wantErr:   true,
			wantErrIs: ErrAccessCheckFailed,
		},
		{
			name:          "manager closed",
			clusterName:   "",
			user:          validUser,
			check:         &AccessCheck{Verb: "get", Resource: "pods"},
			closedManager: true,
			wantErr:       true,
			wantErrIs:     ErrManagerClosed,
		},
		{
			name:        "evaluation error in result",
			clusterName: "",
			user:        validUser,
			check:       &AccessCheck{Verb: "get", Resource: "customresources", APIGroup: "custom.io"},
			setupClient: func(cs *fake.Clientset) {
				cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed:         false,
							EvaluationError: "unable to find resource",
						},
					}, nil
				})
			},
			wantAllowed: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake clients
			scheme := runtime.NewScheme()
			_ = authorizationv1.AddToScheme(scheme)

			fakeClient := fake.NewClientset()
			if tt.setupClient != nil {
				tt.setupClient(fakeClient)
			}

			fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

			provider := &testClientProvider{
				clientset:     fakeClient,
				dynamicClient: fakeDynamic,
				restConfig:    &rest.Config{},
				err:           tt.providerErr,
			}

			manager, err := NewManager(provider)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}

			if tt.closedManager {
				_ = manager.Close()
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := manager.CheckAccess(ctx, tt.clusterName, tt.user, tt.check)

			if tt.wantErr {
				if err == nil {
					t.Error("CheckAccess() expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("CheckAccess() error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				return
			}

			if err != nil {
				t.Errorf("CheckAccess() unexpected error: %v", err)
				return
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("CheckAccess() Allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}

			if result.Denied != tt.wantDenied {
				t.Errorf("CheckAccess() Denied = %v, want %v", result.Denied, tt.wantDenied)
			}

			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("CheckAccess() Reason = %q, want %q", result.Reason, tt.wantReason)
			}
		})
	}
}

func TestManager_CheckAccessAllowed(t *testing.T) {
	validUser := &UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers"},
	}

	tests := []struct {
		name        string
		user        *UserInfo
		check       *AccessCheck
		setupClient func(*fake.Clientset)
		wantErr     bool
		wantErrIs   error
	}{
		{
			name:  "allowed",
			user:  validUser,
			check: &AccessCheck{Verb: "get", Resource: "pods", Namespace: "default"},
			setupClient: func(cs *fake.Clientset) {
				cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: true,
						},
					}, nil
				})
			},
			wantErr: false,
		},
		{
			name:  "denied returns error",
			user:  validUser,
			check: &AccessCheck{Verb: "delete", Resource: "pods", Namespace: "production"},
			setupClient: func(cs *fake.Clientset) {
				cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: false,
							Reason:  "no permission",
						},
					}, nil
				})
			},
			wantErr:   true,
			wantErrIs: ErrAccessDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = authorizationv1.AddToScheme(scheme)

			fakeClient := fake.NewClientset()
			if tt.setupClient != nil {
				tt.setupClient(fakeClient)
			}

			fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

			provider := &testClientProvider{
				clientset:     fakeClient,
				dynamicClient: fakeDynamic,
				restConfig:    &rest.Config{},
			}

			manager, err := NewManager(provider)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = manager.CheckAccessAllowed(ctx, "", tt.user, tt.check)

			if tt.wantErr {
				if err == nil {
					t.Error("CheckAccessAllowed() expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("CheckAccessAllowed() error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				return
			}

			if err != nil {
				t.Errorf("CheckAccessAllowed() unexpected error: %v", err)
			}
		})
	}
}

func TestAccessCheck_SARRequestVerification(t *testing.T) {
	// This test verifies that the SAR request is properly constructed
	user := &UserInfo{
		Email:  "test@example.com",
		Groups: []string{"developers", "testers"},
	}

	check := &AccessCheck{
		Verb:        "delete",
		Resource:    "deployments",
		APIGroup:    "apps",
		Namespace:   "production",
		Name:        "my-deployment",
		Subresource: "scale",
	}

	scheme := runtime.NewScheme()
	_ = authorizationv1.AddToScheme(scheme)

	var capturedAction k8stesting.Action
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		capturedAction = action
		return true, &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})

	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	provider := &testClientProvider{
		clientset:     fakeClient,
		dynamicClient: fakeDynamic,
		restConfig:    &rest.Config{},
	}

	manager, err := NewManager(provider)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = manager.CheckAccess(ctx, "", user, check)
	if err != nil {
		t.Fatalf("CheckAccess() error: %v", err)
	}

	// Verify the request was properly constructed
	if capturedAction == nil {
		t.Fatal("No action was captured")
	}

	createAction, ok := capturedAction.(k8stesting.CreateAction)
	if !ok {
		t.Fatalf("Action was not a CreateAction: %T", capturedAction)
	}

	sar, ok := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
	if !ok {
		t.Fatalf("Created object was not a SelfSubjectAccessReview: %T", createAction.GetObject())
	}

	// Verify all fields were passed correctly
	if sar.Spec.ResourceAttributes == nil {
		t.Fatal("ResourceAttributes was nil")
	}

	ra := sar.Spec.ResourceAttributes
	if ra.Verb != check.Verb {
		t.Errorf("Verb = %q, want %q", ra.Verb, check.Verb)
	}
	if ra.Resource != check.Resource {
		t.Errorf("Resource = %q, want %q", ra.Resource, check.Resource)
	}
	if ra.Group != check.APIGroup {
		t.Errorf("Group = %q, want %q", ra.Group, check.APIGroup)
	}
	if ra.Namespace != check.Namespace {
		t.Errorf("Namespace = %q, want %q", ra.Namespace, check.Namespace)
	}
	if ra.Name != check.Name {
		t.Errorf("Name = %q, want %q", ra.Name, check.Name)
	}
	if ra.Subresource != check.Subresource {
		t.Errorf("Subresource = %q, want %q", ra.Subresource, check.Subresource)
	}
}
