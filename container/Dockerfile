FROM ubuntu:bionic

RUN apt-get update && apt-get install -y wget
ENV GOTRACEBACK=all
ENV GOGC=40
RUN mkdir -p /srv/mr-plotter
ADD assets /srv/mr-plotter/assets
ADD plotter.ini /srv/mr-plotter/
ADD entrypoint.sh /
ADD mr-plotter /bin
ADD hardcodecert /bin
ADD setsessionkeys /bin
RUN chmod a+x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
