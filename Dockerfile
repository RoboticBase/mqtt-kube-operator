FROM alpine:latest
MAINTAINER Nobuyuki Matsui <nobuyuki.matsui@gmail.com>

ARG CONTAINER_BINARY

ENV MQTT_USE_TLS "true"
ENV MQTT_TLS_CA_PATH "/etc/mqtt-kube-operator/certs/DST_Root_CA_X3.pem"
ENV MQTT_USERNAME ""
ENV MQTT_PASSWORD ""
ENV MQTT_HOST "mqtt.example.com"
ENV MQTT_PORT "8883"

COPY ./$CONTAINER_BINARY /opt/mqtt-kube-operator
COPY ./certs/DST_Root_CA_X3.pem /etc/mqtt-kube-operator/certs/DST_Root_CA_X3.pem
WORKDIR /opt
ENTRYPOINT ["/opt/mqtt-kube-operator"]
