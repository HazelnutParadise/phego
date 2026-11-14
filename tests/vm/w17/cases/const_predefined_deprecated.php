<?php
echo E_STRICT, "\n";
echo E_STRICT + 1, "\n";
echo "between\n";
function f()
{
    return E_STRICT;
}
echo f(), f(), "\n";
echo FILE_TEXT, " ", FILE_BINARY, "\n";
$x = E_ALL & ~E_STRICT;
echo $x, "\n";
echo E_STRICT
  . "!\n";
