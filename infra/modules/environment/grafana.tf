resource "grafana_folder" "kosmos" {
  count = var.manage_grafana ? 1 : 0

  title = "Kosmos"
  uid   = "kosmos"
}

resource "grafana_dashboard" "kosmos" {
  count = var.manage_grafana ? 1 : 0

  folder    = grafana_folder.kosmos[0].uid
  overwrite = true
  config_json = jsonencode({
    annotations = {
      list = []
    }
    editable             = true
    fiscalYearStartMonth = 0
    graphTooltip         = 1
    panels = [
      {
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        fieldConfig = {
          defaults = {
            color = {
              mode = "palette-classic"
            }
            unit = "reqps"
          }
          overrides = []
        }
        gridPos = {
          h = 8
          w = 8
          x = 0
          y = 0
        }
        id = 1
        options = {
          legend = {
            calcs       = ["lastNotNull"]
            displayMode = "list"
            placement   = "bottom"
          }
          tooltip = {
            mode = "multi"
            sort = "desc"
          }
        }
        targets = [{
          datasource = {
            type = "loki"
            uid  = var.grafana_logs_datasource_uid
          }
          editorMode   = "code"
          expr         = "sum by (http_response_status_code) (rate({service_name=\"kosmos\"} | http_route != \"\" [$__rate_interval]))"
          legendFormat = "{{http_response_status_code}}"
          queryType    = "range"
          refId        = "A"
        }]
        title = "Request rate by status"
        type  = "timeseries"
      },
      {
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        fieldConfig = {
          defaults = {
            color = {
              mode = "palette-classic"
            }
            unit = "ms"
          }
          overrides = []
        }
        gridPos = {
          h = 8
          w = 8
          x = 8
          y = 0
        }
        id = 2
        options = {
          legend = {
            calcs       = ["lastNotNull"]
            displayMode = "list"
            placement   = "bottom"
          }
          tooltip = {
            mode = "multi"
            sort = "desc"
          }
        }
        targets = [{
          datasource = {
            type = "loki"
            uid  = var.grafana_logs_datasource_uid
          }
          editorMode   = "code"
          expr         = "quantile_over_time(0.95, {service_name=\"kosmos\"} | http_route != \"\" | unwrap duration_ms [$__interval])"
          legendFormat = "p95"
          queryType    = "range"
          refId        = "A"
        }]
        title = "Request latency p95"
        type  = "timeseries"
      },
      {
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        fieldConfig = {
          defaults = {
            color = {
              mode = "thresholds"
            }
            thresholds = {
              mode = "absolute"
              steps = [
                { color = "green", value = null },
                { color = "red", value = 1 },
              ]
            }
          }
          overrides = []
        }
        gridPos = {
          h = 8
          w = 8
          x = 16
          y = 0
        }
        id = 3
        options = {
          colorMode   = "value"
          graphMode   = "area"
          justifyMode = "auto"
          orientation = "auto"
          reduceOptions = {
            calcs  = ["lastNotNull"]
            fields = ""
            values = false
          }
          textMode   = "auto"
          wideLayout = true
        }
        targets = [{
          datasource = {
            type = "loki"
            uid  = var.grafana_logs_datasource_uid
          }
          editorMode = "code"
          expr       = "sum(count_over_time({service_name=\"kosmos\", detected_level=\"error\"}[$__range]))"
          queryType  = "instant"
          refId      = "A"
        }]
        title = "Backend errors"
        type  = "stat"
      },
      {
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        gridPos = {
          h = 10
          w = 12
          x = 0
          y = 8
        }
        id = 4
        options = {
          dedupStrategy      = "none"
          enableLogDetails   = true
          prettifyLogMessage = false
          showCommonLabels   = false
          showLabels         = false
          showTime           = true
          sortOrder          = "Descending"
          wrapLogMessage     = true
        }
        targets = [{
          datasource = {
            type = "loki"
            uid  = var.grafana_logs_datasource_uid
          }
          editorMode = "code"
          expr       = "{service_name=\"kosmos\"} |= \"job\""
          queryType  = "range"
          refId      = "A"
        }]
        title = "Background jobs"
        type  = "logs"
      },
      {
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        gridPos = {
          h = 10
          w = 12
          x = 12
          y = 8
        }
        id = 5
        options = {
          dedupStrategy      = "none"
          enableLogDetails   = true
          prettifyLogMessage = false
          showCommonLabels   = false
          showLabels         = false
          showTime           = true
          sortOrder          = "Descending"
          wrapLogMessage     = true
        }
        targets = [{
          datasource = {
            type = "loki"
            uid  = var.grafana_logs_datasource_uid
          }
          editorMode = "code"
          expr       = "{service_name=\"kosmos\", detected_level=\"error\"}"
          queryType  = "range"
          refId      = "A"
        }]
        title = "Recent backend errors"
        type  = "logs"
      },
      {
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        gridPos = {
          h = 8
          w = 24
          x = 0
          y = 18
        }
        id = 6
        options = {
          dedupStrategy      = "none"
          enableLogDetails   = true
          prettifyLogMessage = false
          showCommonLabels   = false
          showLabels         = false
          showTime           = true
          sortOrder          = "Descending"
          wrapLogMessage     = true
        }
        targets = [{
          datasource = {
            type = "loki"
            uid  = var.grafana_logs_datasource_uid
          }
          editorMode = "code"
          expr       = "{app_id=\"902\"} | detected_level=\"error\""
          queryType  = "range"
          refId      = "A"
        }]
        title = "Frontend errors"
        type  = "logs"
      },
    ]
    refresh       = "1m"
    schemaVersion = 41
    tags          = ["kosmos", var.environment, "managed-by-opentofu"]
    templating = {
      list = []
    }
    time = {
      from = "now-6h"
      to   = "now"
    }
    timezone = "browser"
    title    = "Kosmos ${title(var.environment)} Overview"
    uid      = "kosmos-${var.environment}"
    version  = 0
  })
}

resource "grafana_rule_group" "kosmos" {
  count = var.manage_grafana ? 1 : 0

  name             = "Kosmos ${title(var.environment)}"
  folder_uid       = grafana_folder.kosmos[0].uid
  interval_seconds = 60

  rule {
    name           = "Kosmos frontend error detected"
    for            = "0s"
    condition      = "B"
    no_data_state  = "OK"
    exec_err_state = "Alerting"
    is_paused      = false

    annotations = {
      description = "Faro recorded a production browser error. Check the frontend error panel and correlated trace in the Kosmos dashboard."
      summary     = "Kosmos frontend error detected"
    }

    labels = {
      environment = var.environment
      service     = "kosmos"
      severity    = "error"
    }

    data {
      ref_id         = "A"
      query_type     = "range"
      datasource_uid = var.grafana_logs_datasource_uid

      relative_time_range {
        from = 300
        to   = 0
      }

      model = jsonencode({
        datasource = {
          type = "loki"
          uid  = var.grafana_logs_datasource_uid
        }
        editorMode    = "code"
        expr          = "sum(count_over_time({app_id=\"902\", detected_level=\"error\"}[5m]))"
        intervalMs    = 1000
        maxDataPoints = 43200
        queryType     = "range"
        refId         = "A"
      })
    }

    data {
      ref_id         = "B"
      datasource_uid = "-100"

      relative_time_range {
        from = 0
        to   = 0
      }

      model = jsonencode({
        conditions = [{
          evaluator = {
            params = [0]
            type   = "gt"
          }
          operator = {
            type = "and"
          }
          query = {
            params = ["A"]
          }
          reducer = {
            params = []
            type   = "last"
          }
          type = "query"
        }]
        datasource = {
          type = "__expr__"
          uid  = "-100"
        }
        hide          = false
        intervalMs    = 1000
        maxDataPoints = 43200
        refId         = "B"
        type          = "classic_conditions"
      })
    }
  }
}
