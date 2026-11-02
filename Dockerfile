# Build the SPA, embed it into a static Go binary, run it as non-root.

FROM node:24.22.2-alpine3.24 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26.10-alpine3.24 AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/internal/spa/dist ./internal/spa/dist
RUN go build -trimpath -ldflags="-s -w" -o /out/moimroom ./cmd/moimroom

# distroless static: CA certificates, no shell, uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/moimroom /moimroom
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/moimroom"]
