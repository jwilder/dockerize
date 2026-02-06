FROM ubuntu:noble

SHELL ["/bin/bash", "-Eeuxo", "pipefail", "-c"]

RUN <<EOF
    apt-get update && apt-get install -y wget nginx

    echo "daemon off;" >> /etc/nginx/nginx.conf
    ln -sf /dev/stdout /var/log/nginx/access.log
    ln -sf /dev/stderr /var/log/nginx/error.log
EOF

RUN wget https://github.com/invarnt/dockerize/releases/download/v0.1.0/dockerize-linux-amd64-v0.1.0.tar.gz \
    && tar -C /usr/local/bin -xvzf dockerize-linux-amd64-v0.1.0.tar.gz

ADD default.tmpl /etc/nginx/sites-available/default.tmpl

EXPOSE 80

CMD ["dockerize", "-template", "/etc/nginx/sites-available/default.tmpl:/etc/nginx/sites-available/default", "nginx"]
