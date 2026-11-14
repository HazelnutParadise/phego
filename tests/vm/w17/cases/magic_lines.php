<?php
echo
  __LINE__,
  "\n";
$a = [
    __LINE__,
    __LINE__
];
echo $a[0], $a[1], "\n";
echo __LINE__ .
  __LINE__ .
  __LINE__, "\n";
function f(
    $l = __LINE__
) {
    return $l . __LINE__;
}
echo f(), "\n";
