# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang:latest

# Copy the local package files to the container's workspace.
ADD . /go/src/github.com/agile-leaf/50mm

# get the binary together
WORKDIR /go/src/github.com/agile-leaf/50mm
RUN go get -v


# get the deploy folder structure in working condition
RUN mkdir /deploy
WORKDIR /deploy

# add in the files we need in the workdir
RUN mv /go/bin/50mm .
ADD static ./static
ADD templates ./templates
RUN mkdir config

# get all the working parts in place to get running
ENV FIFTYMM_PORT=80
ENV FIFTYMM_CONFIG_DIR=/deploy/config

# Run the outyet command by default when the container starts.
CMD /deploy/50mm

# Document that the service listens on port 80.
EXPOSE 80