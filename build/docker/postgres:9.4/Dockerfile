FROM postgres:9.4

COPY --chown=postgres postgresql.conf /usr/share/postgresql/

# Check with $ psql -U mmuser mattermost_test -c 'SHOW config_file'
CMD ["postgres", "-c", "config_file=/usr/share/postgresql/postgresql.conf"]
