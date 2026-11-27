<?php
$e = new
  Exception(
    "m"
  );
echo $e->getLine(), "\n";
$f = new Exception(
  "a" .
  "b"
);
echo $f->getLine(), "\n";
function g() {
  return new Exception(
    "z");
}
echo g()->getLine(), " ", g()->getTrace()[0]["line"], "\n";
