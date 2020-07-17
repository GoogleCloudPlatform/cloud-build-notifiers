# Cloud Build BigQuery Notifier

This notifier pushes build data to a BigQuery instance.

*Alpha Feature - Under Development*

SQL queries through the BigQuery CLI:

bq query '\<SQL QUERY\>'

Legacy SQL dialect is set on default for the BigQuery CLI and must be disabled for the example queries to work.
This can be done by editing ~/.bigqueryrc and adding the following lines:

```
[query]
--use_legacy_sql=false
```

Example Queries:
```
# Listing overall build history

SELECT * FROM `dataset.table`

# Aggregating Build Counts by Status

SELECT STATUS,  COUNT(*) FROM `dataset.table` GROUP BY STATUS

# Getting Daily Deployment Frequency for Current Week

SELECT DAY, COUNT(STATUS) AS Deployments FROM (SELECT DATETIME_TRUNC(CreateTIme, WEEK) AS WEEK, DATETIME_TRUNC(CreateTime, DAY) AS DAY, STATUS FROM `dataset.table` WHERE STATUS="SUCCESS") WHERE WEEK = DATETIME_TRUNC(CURRENT_DATETIME(), WEEK) GROUP BY DAY'

# Calculating BuildTimes

SELECT CreateTime, DATETIME_DIFF(FinishTime, StartTime, SECOND) as BuildTime FROM `dataset.table`  WHERE STATUS = "SUCCESS" ORDER BY BuildTime

# Getting Build Statuses for the Current Day

SELECT DAY, STATUS FROM (SELECT DATETIME_TRUNC(CreateTime, DAY) AS DAY, STATUS FROM `dataset.table`) WHERE DAY = DATETIME_TRUNC(CURRENT_DATETIME(), DAY)
```
