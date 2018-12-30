FROM ubuntu:18.04
MAINTAINER Jason Wilder mail@jasonwilder.com

# Install Nginx.
RUN apt-get update && apt-get install -y wget nginx

RUN wget https://github.com/jwilder/dockerize/releases/download/v0.6.1/dockerize-linux-amd64-v0.6.1.tar.gz \
    && tar -C /usr/local/bin -xvzf dockerize-linux-amd64-v0.6.1.tar.gz

RUN echo "daemon off;" >> /etc/nginx/nginx.conf

ADD default.tmpl /etc/nginx/sites-available/default.tmpl

EXPOSE 80

CMD ["dockerize", "-template", "/etc/nginx/sites-available/default.tmpl:/etc/nginx/sites-available/default", "-stdout", "/var/log/nginx/access.log", "-stderr", "/var/log/nginx/error.log", "nginx"]
