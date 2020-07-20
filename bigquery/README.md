# Cloud Build BigQuery Notifier

This notifier pushes build data to a BigQuery instance.

*Alpha Feature - Under Development*

## Accessing build insights with SQL queries through the BigQuery CLI:

To access BQ data through queries, run the following command below.


```bash
$ bq query '<SQL QUERY>'
```
More detailed information can be found here: [BQ CLI Reference](https://cloud.google.com/bigquery/docs/bq-command-line-tool)

Legacy SQL dialect is set on default for the BigQuery CLI and must be disabled for the example queries to work.
This can be done by adding the the ```--nouse_legacy_sql``` flag:

```bash
$ bq query --nouse_legacy_sql '<SQL QUERY>'
```

Alternatively, removing the flag requirement would require editing ```~/.bigqueryrc``` and adding the following lines:

```
[query]
--use_legacy_sql=false
```
More information can be found here: [Switching SQL Dialects](https://cloud.google.com/bigquery/docs/reference/standard-sql/enabling-standard-sql).

### Example Queries:

```sql
# Listing overall build history

SELECT * FROM `dataset.table`

# Aggregating build counts by status

SELECT STATUS, COUNT(*) 
FROM `dataset.table` 
GROUP BY STATUS

# Getting daily deployment frequency for current week

SELECT DAY, COUNT(STATUS) AS Deployments 
FROM (SELECT DATETIME_TRUNC(CreateTime, WEEK) AS WEEK, 
      DATETIME_TRUNC(CreateTime, DAY) AS DAY, 
      STATUS 
      FROM `dataset.table` 
      WHERE STATUS="SUCCESS") 
WHERE WEEK = DATETIME_TRUNC(CURRENT_DATETIME(), WEEK) 
GROUP BY DAY

# Calculating build times

SELECT CreateTime, DATETIME_DIFF(FinishTime, StartTime, SECOND) as BuildTime 
FROM `dataset.table`  
WHERE STATUS = "SUCCESS" 
ORDER BY BuildTime

# Getting build statuses for the current day

SELECT DAY, STATUS 
FROM (SELECT DATETIME_TRUNC(CreateTime, DAY) AS DAY, 
      STATUS FROM `dataset.table`) 
WHERE DAY = DATETIME_TRUNC(CURRENT_DATETIME(), DAY)
```
