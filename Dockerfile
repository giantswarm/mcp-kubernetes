FROM gsoci.azurecr.io/giantswarm/alpine:3.20.3-giantswarm
FROM scratch

COPY --from=0 /etc/passwd /etc/passwd
COPY --from=0 /etc/group /etc/group

ADD mcp-kubernetes /
USER giantswarm

ENTRYPOINT ["/mcp-kubernetes"]
