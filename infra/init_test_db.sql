-- init-test-db.sql
CREATE USER testuser WITH PASSWORD 'testpass';
CREATE DATABASE testdb;
GRANT ALL PRIVILEGES ON DATABASE testdb TO testuser;
ALTER DATABASE testdb OWNER TO testuser;