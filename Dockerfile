# Use a minimal base image
FROM alpine:latest

# Create a file with dummy secret keys
RUN echo "secret_key=ASIAJ73N6GYZRLJCM52Q" > /dummy_secrets.txt && \
    echo "api_token=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A" >> /dummy_secrets.txt

RUN echo "ASIAIQAP7NCOV4IOP6HQ"
# Print the contents of the file during the build
RUN cat /dummy_secrets.txt

RUN cp /dummy_secres.txt . 