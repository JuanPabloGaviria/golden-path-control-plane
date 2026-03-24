FROM golang:1.25 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG APP_BIN=api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./cmd/${APP_BIN}

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/app /app

USER nonroot:nonroot

ENTRYPOINT ["/app"]
