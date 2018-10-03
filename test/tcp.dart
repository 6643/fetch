import 'dart:io';
import 'dart:convert';

main(List<String> args) {
  // _get();
  _post();
}

_get() {
  Socket.connect('httpbin.org', 80).then((tcp) {
    print(tcp);
    tcp.listen((onData) {
      print(utf8.decode(onData));
    });
    tcp.write(
        'GET /get HTTP/1.1\r\nHost: httpbin.org\r\naccept: application/json\r\nConnection: close\r\n\r\n');
  }).catchError(() {
    print('onError');
  });
}

_post() {
  Socket.connect('httpbin.org', 80).then((tcp) {
    print(tcp);
    tcp.listen((onData) {
      print(utf8.decode(onData));
      tcp.close();
    });

    tcp.write('POST /post?a=b&a=c HTTP/1.1\r\n');
    tcp.write('Host: httpbin.org\r\n');
    tcp.write('Content-Type: application/json;charset=utf-8\r\n');
    tcp.write('Content-Length: 9\r\n');
    tcp.write('Connection: close\r\n\r\n');
    tcp.write('{"a":"b"}');

    tcp.flush();
  }).catchError(() {
    print('onError');
  });
}
