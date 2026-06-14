# Slim since we don't need extra build tools and Alpine might break clip
FROM python:3.12-slim

WORKDIR /app

ENV PYTHONUNBUFFERED=1

# Install dependencies first for layer caching.
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Copy the source.
COPY src/ ./src/

ENV PYTHONPATH=/app/src

CMD ["python", "-m", "worker.main"]