FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH
COPY dist/kosmos_linux_${TARGETARCH}*/kosmos /kosmos
COPY frontend/dist /web
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/kosmos"]
