FROM nginx:1.9
MAINTAINER Jason Wilder mail@jasonwilder.com

# RUN wget https://github.com/markriggins/dockerize/releases/download/v0.2.0/dockerize-linux-amd64-v0.0.4.tar.gz
# RUN tar -C /usr/local/bin -xvzf dockerize-linux-amd64-v0.2.0.tar.gz

COPY .dockerize_linux_amd64.tar.gz /tmp/
RUN  cd /usr/local/bin && \
    tar -xzv --strip-components=1 -f /tmp/.d*.gz dockerize_linux_amd64/dockerize && \
    rm /tmp/.d*.gz

COPY default.tmpl /etc/nginx/sites-available/default.tmpl
COPY overlays /tmp/overlays

EXPOSE 80

ENV DEPLOYMENT_ENV=staging

#  These options do not work well on virtualbox osx -- it pegs one CPU
#     "-poll", \
#     "-stdout", "/var/log/nginx/access.log", \
#     "-stderr", "/var/log/nginx/error.log", \

CMD [ "dockerize", \
     "-template", "/etc/nginx/sites-available/default.tmpl:/etc/nginx/sites-available/default", \
     "-overlay", "/tmp/overlays/_common/html:/usr/share/nginx/", \
     "-overlay", "/tmp/overlays/{{ .Env.DEPLOYMENT_ENV }}/html:/usr/share/nginx/", \
     "-poll", \
     "-stdout", "/var/log/nginx/access.log", \
     "-stderr", "/var/log/nginx/error.log", \
     "--", "nginx", "-g", "daemon off;"]
