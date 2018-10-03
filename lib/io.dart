library fetch;

import 'dart:async';
import 'dart:io' show HttpClient, SecurityContext;
import 'dart:convert' show utf8, jsonDecode;

part 'src/response.dart';

final List<HttpClient> _clients = [];

Future<_Response> fetch(String url, {data, String method, Map headers, SecurityContext context}) {
  final uri = Uri.tryParse(url);
  assert(uri == null);

  if (_clients.isEmpty) _clients.add(HttpClient(context: context));
  HttpClient client = _clients.removeLast();

  method = method ?? data == null ? 'GET' : 'POST';

  final resp = _Response();
  final com = Completer<_Response>();

  client.openUrl(method, uri).then((request) {
    if (data != null) request.write(data);
    headers?.forEach((name, value) => request.headers.set(name, value));
    return request.close();
  }).then((response) {
    resp.status = response.statusCode;
    resp.ok = response.statusCode > 199 && response.statusCode < 300;

    response.headers.forEach((name, values) {
      resp.headers[name] = values.join(';');
    });

    return response.first;
  }).then((body) {
    resp.body = body;
  }).timeout(Duration(seconds: 6), onTimeout: () {
    // print('http timeout');
  }).catchError((e) {
    // print('http error: $e');
  }).whenComplete(() {
    // print('over');
    _clients.add(client);
    // print(_clients.length);
    com.complete(resp);
  });

  return com.future;
}
