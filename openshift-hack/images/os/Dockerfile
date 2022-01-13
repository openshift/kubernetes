FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.17-openshift-4.10 AS build

COPY --from=registry.ci.openshift.org/ocp/4.10:machine-os-content /srv/ /srv/
COPY --from=registry.ci.openshift.org/ocp/4.10:machine-os-content /extensions/ /extensions/
WORKDIR /
COPY install.sh .
RUN ./install.sh

FROM scratch
COPY --from=build /srv/ /srv/
COPY --from=build /extensions/ /extensions/

LABEL io.openshift.build.version-display-names="machine-os=rhcos image for testing openshift kubernetes kubelet only- if you see this outside of PR runs for openshift kubernetes- you found an urgent blocker bug" \
      io.openshift.build.versions="machine-os=1.2.3-testing-if-you-see-this-outside-of-PR-runs-for-openshift-kubernetes-you-found-an-urgent-blocker-bug"
