FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o relay .

FROM alpine:3.19
COPY --from=build /app/relay /relay
CMD ["/relay"]
