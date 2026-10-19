FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nvuto-consent ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nvuto-consent /nvuto-consent
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/nvuto-consent"]
