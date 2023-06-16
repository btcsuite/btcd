name: Docker images build and publish

on:
  push:
    tags:
      - v*
  # Allows you to run this workflow manually from the Actions tab
  workflow_dispatch:

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}
  # Build for default OS, linux, and common CPU architectures
  # Reference https://github.com/docker/setup-buildx-action#quick-start
  TPLATFORMS: linux/amd64,linux/arm64,linux/arm,linux/386

jobs:
  build-push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout repository
        uses: actions/checkout@v2

      - name: Docker Setup Buildx
        id: buildx
        uses: docker/setup-buildx-action@94ab11c41e45d028884a99163086648e898eed25

      - name: Log in to the Container registry
        uses: docker/login-action@f054a8b539a109f9f41c372932f1ae047eff08c9
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata (tags, labels) for Docker
        id: meta
        uses: docker/metadata-action@98669ae865ea3cffbcbaa878cf57c20bbf1c6c38
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
       
      - name: Build and push Docker images
        uses: docker/build-push-action@ac9327eae2b366085ac7f6a2d02df8aa8ead720a
        with:
          file: .github/workflows/Dockerfile
          labels: ${{ steps.meta.outputs.labels }}
          platforms: ${{ env.TPLATFORMS }}
          push: true
          tags: ${{ steps.meta.outputs.tags }}
