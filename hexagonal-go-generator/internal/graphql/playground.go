// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// playground.go - serves a single-page GraphiQL UI bound to /graphql.
package graphql

import "net/http"

// PlaygroundHandler serves GraphiQL.
func PlaygroundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(graphiqlHTML))
	}
}

const graphiqlHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Hexagonal Go - GraphiQL</title>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3.0.10/graphiql.min.css">
  <style>body { margin: 0; }</style>
</head>
<body>
  <div id="graphiql" style="height: 100vh;"></div>
  <script src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script src="https://unpkg.com/graphiql@3.0.10/graphiql.min.js"></script>
  <script>
    ReactDOM.render(
      React.createElement(GraphiQL, { fetcher: (params) =>
        fetch('/graphql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(params),
        }).then(r => r.json())
      }),
      document.getElementById('graphiql'),
    );
  </script>
</body>
</html>`
