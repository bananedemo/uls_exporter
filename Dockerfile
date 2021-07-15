FROM golang
WORKDIR /build
COPY . .
RUN go build -v .

FROM gcr.io/distroless/base
COPY --from=0 /build/uls_exporter /bin/uls_exporter
CMD ["/bin/uls_exporter"]