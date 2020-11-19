# Cloud Build BigQuery Notifier

This notifier pushes build data to a BigQuery instance.

## Dataset and table setup

The BQ notifier initializes datasets and tables automatically.
Dataset identifiers in the BigQuery Notifier config can refer to existing or nonexistent datasets. 

Table identifiers in the BigQuery Notifier config can refer to either:
1. A nonexistent table (will be created upon deployment of the notifier)
2. An empty table not yet initialized with a schema.
3. An existing table with a schema that matches the bq notifier schema specifications.

References to already existing tables with differing schemas will throw errors upon writing.

## Accessing build insights with SQL queries through the BigQuery CLI:

To access BQ data through queries, run the following command below.

```bash
$ bq query '<SQL QUERY>'
```
Adding the `--format=prettyjson` flag allows for more readable output.

More detailed information can be found here: [BQ CLI Reference](https://cloud.google.com/bigquery/docs/bq-command-line-tool)

Legacy SQL dialect is set on default for the BigQuery CLI and must be disabled for the example queries to work.
This can be done by adding the the `--nouse_legacy_sql` flag:

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

SELECT * FROM `projectID.datasetName.tableName`

# Aggregating build counts by status

SELECT STATUS, COUNT(*) 
FROM `projectID.datasetName.tableName`
GROUP BY STATUS

# Getting daily deployment frequency for current week

SELECT DAY, COUNT(STATUS) AS Deployments 
FROM (SELECT DATETIME_TRUNC(CreateTime, WEEK) AS WEEK, 
      DATETIME_TRUNC(CreateTime, DAY) AS DAY, 
      STATUS 
      FROM `projectID.datasetName.tableName` 
      WHERE STATUS="SUCCESS") 
WHERE WEEK = DATETIME_TRUNC(CURRENT_DATETIME(), WEEK) 
GROUP BY DAY

# Calculating build times

SELECT CreateTime, DATETIME_DIFF(FinishTime, StartTime, SECOND) as BuildTime 
FROM `projectID.datasetName.tableName`  
WHERE STATUS = "SUCCESS" 
ORDER BY BuildTime

# Getting build statuses for the current day

SELECT DAY, STATUS 
FROM (SELECT DATETIME_TRUNC(CreateTime, DAY) AS DAY, 
      STATUS FROM `projectID.datasetName.tableName`) 
WHERE DAY = DATETIME_TRUNC(CURRENT_DATETIME(), DAY)
```
