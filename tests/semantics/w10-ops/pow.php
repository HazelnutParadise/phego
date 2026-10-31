<?php
// Reads "x y" pairs of 16-hex-digit IEEE-754 bit patterns, one per line, and
// prints the bits of x ** y as computed by PHP.
set_error_handler(fn() => true); // deprecations do not change the result
while (($line = fgets(STDIN)) !== false) {
    [$x, $y] = explode(' ', rtrim($line, "\n"));
    $r = unpack('E', hex2bin($x))[1] ** unpack('E', hex2bin($y))[1];
    echo bin2hex(pack('E', $r)), "\n";
}
