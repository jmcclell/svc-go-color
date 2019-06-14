docker build -t colro-dev -f docker/Dockerfile-dev
ocker run --rm -it -p 8000:3000 -v $(PWD):/go/src/app --network dev color-dev

