.PHONY : build run-staging run-prod

build: 
	cd ../..; goxc -os linux -arch amd64
	cp -f $(shell bash -xc 'find $(GOPATH)/bin -name dockerize_linux_amd64.tar.gz| head -1') .dockerize_linux_amd64.tar.gz
	docker build -t dockerized-nginx .
	rm .dockerize_linux_amd64.tar.gz


run-staging: 
	docker rm -f dockerized-nginx 2>/dev/null || true
	docker run -d -p 80:80 -e DEPLOYMENT_ENV=staging --name dockerized-nginx dockerized-nginx
	open http://$(shell docker-machine ip $(shell docker-machine active))/robots.txt

run-prod: 
	docker rm -f dockerized-nginx 2>/dev/null || true
	docker run -d -p 80:80 -e DEPLOYMENT_ENV=prod --name dockerized-nginx dockerized-nginx
	open http://$(shell docker-machine ip $(shell docker-machine active))/robots.txt
