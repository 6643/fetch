class Request {
  String method;
  String url;
  dynamic data;

  Map headers;
  Request(this.method, this.url, this.data, this.headers);
}
