# Build the Go service. Java 21 is not needed: the service is Go (net/http),
# a deliberate, documented deviation from the Spring Boot spec. See REPORT.md.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /service .

FROM gcr.io/distroless/static-debian12
COPY --from=build /service /service
EXPOSE 8443
ENTRYPOINT ["/service"]
