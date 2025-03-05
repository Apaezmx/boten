# Stage 1: Builder stage
FROM --platform=linux/amd64 golang:1.22-alpine
WORKDIR /app
COPY . ./
RUN go mod download
RUN go build -o myapp
ENV GOOGLE_APPLICATION_CREDENTIALS=/app/retaissance-7dfc39aaeecc.json
EXPOSE 8080
CMD ["./myapp"]