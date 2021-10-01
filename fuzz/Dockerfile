FROM golang:1.16-buster as builder

RUN apt-get update && apt-get install -y clang git
RUN git clone --branch fuzz1 --depth 1 https://github.com/AdamKorcz/helm-controller /helm-controller
COPY fuzz.go /helm-controller/controllers/
RUN cd /helm-controller/controllers \
	&& go get github.com/AdaLogics/go-fuzz-headers

RUN cd / \
	&& go get -u github.com/mdempsky/go114-fuzz-build

RUN mkdir /fuzzers
RUN cd /helm-controller/controllers \
	&& go114-fuzz-build -o FuzzHelmreleaseComposeValues.a -func FuzzHelmreleaseComposeValues . \
	&& clang -o /fuzzers/FuzzHelmreleaseComposeValues FuzzHelmreleaseComposeValues.a -fsanitize=fuzzer
RUN cd /helm-controller/controllers \
	&& go114-fuzz-build -o FuzzHelmreleasereconcile.a -func FuzzHelmreleasereconcile . \
	&& clang -o /fuzzers/FuzzHelmreleasereconcile FuzzHelmreleasereconcile.a -fsanitize=fuzzer

# Uncomment here to run the fuzzer
#RUN /fuzzers/FuzzHelmreleaseComposeValues
