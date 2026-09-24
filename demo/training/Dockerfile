FROM python:3.13-slim
WORKDIR /app
COPY train.py /app/train.py
USER 65534:65534
ENTRYPOINT ["python", "-u", "/app/train.py"]
