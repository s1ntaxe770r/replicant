FROM golang:latest as builder

WORKDIR /app

COPY . . 

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server main.go

FROM alpine 

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/server /server

EXPOSE 8080 

ENTRYPOINT ["/server"]
