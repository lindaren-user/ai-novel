FROM golang:1.25-alpine AS builder

WORKDIR /app

# 国内需要
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off 

COPY be/go.mod be/go.sum ./
RUN go mod download

COPY be/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/ai-novel-server ./cmd/server

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/ai-novel-server /app/ai-novel-server
COPY be/internal/service/ai_skills /app/ai_skills

EXPOSE 8080

CMD ["/app/ai-novel-server"]
