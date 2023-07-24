FROM quay.io/astronomer/:new-version
USER root
RUN pip install pip-tools
RUN pip freeze > req.txt
RUN cat requirements.txt >> req.txt
RUN sed -i '/\.whl/d' req.txt
RUN python -m piptools compile --verbose req.txt -o conflict-test-results.txt
