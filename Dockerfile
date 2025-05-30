FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./main.go

FROM alpine:latest
RUN apk update
RUN apk add blender curl py3-pip
RUN pip install --root-user-action ignore --break-system-packages numpy scipy trimesh
RUN apk del py3-pip

WORKDIR /root/
COPY --from=builder /app/server .
COPY main.py.tmpl .
EXPOSE 1212
CMD ["./server"]
