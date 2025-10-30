# ========= GO =========
FROM golang:1.25-alpine AS buildergo

WORKDIR /app
COPY ./ ./
RUN go build -o /app/helheim-master ./cmd/helheim-master/main.go
RUN go build -o /app/helheim-slave ./cmd/helheim-slave/main.go


# ========= APP =========
FROM ubuntu:22.04

# timezone
ARG TZ=UTC
ENV TZ ${TZ}
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# user
ARG PUID=1000
ENV PUID ${PUID}
ARG PGID=1000
ENV PGID ${PGID}

RUN groupadd -g $PGID app && \
  useradd app -u $PUID -g $PGID -m -s /bin/bash

# files
WORKDIR /app
COPY --from=buildergo /app/helheim-master /app/helheim-master
COPY --from=buildergo /app/helheim-slave /app/helheim-slave
RUN mkdir -p /var/sync && chown -R $PUID:$PGID /var/sync

# run
USER app