FROM golang:1.17 as builder

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/ydb-kubernetes-operator/main.go

FROM alpine:3.15
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
