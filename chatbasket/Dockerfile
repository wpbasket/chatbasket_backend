FROM golang:1.24.4-alpine AS build_base
RUN apk add --no-cache git
WORKDIR /app

COPY . .
RUN go mod download

RUN GOOS=linux CGO_ENABLED=0 go build -o bootstrap ./app
FROM alpine:3.22.0
RUN apk add ca-certificates

# To use the AWS Lambda Web Adapter in the docker image
COPY --from=public.ecr.aws/awsguru/aws-lambda-adapter:0.9.1 /lambda-adapter /opt/extensions/lambda-adapter
COPY --from=build_base /app/bootstrap /app/bootstrap

ENV PORT=8000 AWS_LWA_ASYNC_INIT=true
EXPOSE 8000

CMD ["/app/bootstrap"]