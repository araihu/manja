package render

type RequestSampleTarget struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Target   string `json:"target"`
	Client   string `json:"client"`
	Language string `json:"language"`
}

func RequestSampleTargets() []RequestSampleTarget {
	return []RequestSampleTarget{
		{Value: "shell:curl", Label: "Shell / cURL", Target: "shell", Client: "curl", Language: "shell"},
		{Value: "shell:httpie", Label: "Shell / HTTPie", Target: "shell", Client: "httpie", Language: "shell"},
		{Value: "shell:wget", Label: "Shell / Wget", Target: "shell", Client: "wget", Language: "shell"},
		{Value: "javascript:fetch", Label: "JavaScript / fetch", Target: "javascript", Client: "fetch", Language: "javascript"},
		{Value: "javascript:axios", Label: "JavaScript / Axios", Target: "javascript", Client: "axios", Language: "javascript"},
		{Value: "node:fetch", Label: "Node.js / fetch", Target: "node", Client: "fetch", Language: "javascript"},
		{Value: "python:requests", Label: "Python / Requests", Target: "python", Client: "requests", Language: "python"},
		{Value: "go:native", Label: "Go / NewRequest", Target: "go", Client: "native", Language: "go"},
		{Value: "java:okhttp", Label: "Java / OkHttp", Target: "java", Client: "okhttp", Language: "java"},
		{Value: "php:curl", Label: "PHP / cURL", Target: "php", Client: "curl", Language: "php"},
		{Value: "ruby:native", Label: "Ruby / net::http", Target: "ruby", Client: "native", Language: "ruby"},
		{Value: "csharp:restsharp", Label: "C# / RestSharp", Target: "csharp", Client: "restsharp", Language: "csharp"},
		{Value: "powershell:webrequest", Label: "Powershell / Invoke-WebRequest", Target: "powershell", Client: "webrequest", Language: "powershell"},
		{Value: "powershell:restmethod", Label: "Powershell / Invoke-RestMethod", Target: "powershell", Client: "restmethod", Language: "powershell"},
		{Value: "swift:urlsession", Label: "Swift / URLSession", Target: "swift", Client: "urlsession", Language: "swift"},
		{Value: "kotlin:okhttp", Label: "Kotlin / OkHttp", Target: "kotlin", Client: "okhttp", Language: "kotlin"},
		{Value: "http:http1.1", Label: "HTTP / HTTP/1.1", Target: "http", Client: "http1.1", Language: "http"},
		{Value: "c:libcurl", Label: "C / Libcurl", Target: "c", Client: "libcurl", Language: "c"},
		{Value: "clojure:clj_http", Label: "Clojure / clj-http", Target: "clojure", Client: "clj_http", Language: "clojure"},
		{Value: "objc:nsurlsession", Label: "Objective-C / NSURLSession", Target: "objc", Client: "nsurlsession", Language: "objc"},
		{Value: "ocaml:cohttp", Label: "OCaml / CoHTTP", Target: "ocaml", Client: "cohttp", Language: "ocaml"},
		{Value: "r:httr", Label: "R / httr", Target: "r", Client: "httr", Language: "r"},
	}
}
