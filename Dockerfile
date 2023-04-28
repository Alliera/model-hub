FROM golang:1.19.1-bullseye

WORKDIR /src

# download dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# build and install clerk executable
COPY . .
RUN go build -ldflags "-s -w" -o /model-hub

FROM huggingface/transformers-pytorch-gpu:latest

WORKDIR /bin
COPY --from=0 /model-hub /bin/model-hub
COPY ./worker.py /bin/worker.py

CMD model-hub
