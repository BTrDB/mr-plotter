FROM ubuntu:xenial


RUN apt-get update && apt-get install -y  wget git
ENV GOTRACEBACK=all
ENV GOGC=40
RUN cd /srv && \
    git clone https://github.com/BTrDB/mr-plotter.git && \
    cd mr-plotter
ADD entrypoint.sh /
ADD mr-plotter /bin
ADD hardcodecert /bin
ADD setsessionkeys /bin
RUN chmod a+x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
