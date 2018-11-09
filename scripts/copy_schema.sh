#!/bin/sh

set -xe

mv ../docker/sql/schema.sql ../docker/sql/schema.old.sql
cp ../db/schema.sql ../docker/sql/schema.sql
