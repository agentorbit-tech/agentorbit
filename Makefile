.PHONY: web build-processing build-proxy

web:
	cd web && npm install && npm run build
	rm -rf processing/web/dist
	mkdir -p processing/web/dist
	cp -r web/dist/. processing/web/dist/
	touch processing/web/dist/.gitkeep

build-processing: web
	cd processing && go build -o ../bin/processing ./cmd/processing

build-proxy:
	cd proxy && go build -o ../bin/proxy ./cmd/proxy
