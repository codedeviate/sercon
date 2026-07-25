// text.case — convert identifiers between naming conventions.
const id = "getHTTPResponseCode";

runtime.assert.equal(text.case.snake(id), "get_http_response_code");
runtime.assert.equal(text.case.kebab(id), "get-http-response-code");
runtime.assert.equal(text.case.pascal(id), "GetHttpResponseCode");
runtime.assert.equal(text.case.camel("get-http-response-code"), "getHttpResponseCode");
runtime.assert.equal(text.case.screamingSnake(id), "GET_HTTP_RESPONSE_CODE");
runtime.assert.equal(text.case.dot(id), "get.http.response.code");

// Dynamic dispatch + the primitives.
runtime.assert.equal(text.case.convert(id, "title"), "Get Http Response Code");
runtime.assert.equal(JSON.stringify(text.case.split(id)), JSON.stringify(["get","http","response","code"]));
runtime.assert.equal(text.case.detect("get_http"), "snake");

runtime.log("text.case:", text.case.names().length, "cases");
for (const name of text.case.names()) {
  runtime.log(`  ${name.padEnd(16)} ${text.case.convert(id, name)}`);
}
