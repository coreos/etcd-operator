FROM golang:1.9

RUN curl -L https://github.com/golang/dep/releases/download/v0.3.2/dep-linux-amd64 -o /usr/local/bin/dep \
    && chmod +x /usr/local/bin/dep \
    && go get honnef.co/go/tools/cmd/gosimple \
    && go get honnef.co/go/tools/cmd/unused
