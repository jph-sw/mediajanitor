# goreleaser passes a pre-built binary into this build context.
# This stage exists only to supply ca-certificates to the scratch image.
FROM alpine:3.19 AS certs
RUN apk --no-cache add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY mediajanitor /mediajanitor
ENTRYPOINT ["/mediajanitor"]
