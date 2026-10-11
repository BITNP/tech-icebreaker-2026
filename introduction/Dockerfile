# 两段构建：node 出前端静态产物，go 把产物嵌进单个二进制。
FROM node:22-alpine AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY babel.config.js jsconfig.json vue.config.js ./
COPY src ./src
COPY public ./public
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/dist ./dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/icebreaker ./cmd/icebreaker

FROM alpine
WORKDIR /app
COPY --from=build /out/icebreaker /app/icebreaker
# config.yaml 与 data/*.db 都不进镜像，部署时从宿主机挂载到 /app/config.yaml 和 /app/data。
RUN mkdir -p /app/data
EXPOSE 8080
ENTRYPOINT ["/app/icebreaker", "-config", "/app/config.yaml"]
