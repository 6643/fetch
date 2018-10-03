part of fetch;

class _Response {
  List<int> body;
  Map headers = {};
  bool ok = false;
  int status = 0;

  _Response();

  Map args() {}
  Map form() {}
  Map files() {}
  dynamic json() => jsonDecode(text());
  String text() => utf8.decode(body);
}
