<?php
// CVE: CVE-2017-6441
// A crafted declare(ticks=...) value reached _zval_get_long_func_ex as a
// NULL value. PHP 8.5 takes any literal and converts it to an integer,
// warning for a float outside the integer range. The cases ending in
// _null, _true and _constant cover the forms it rejects.
declare(ticks="1");
declare(ticks="abc");
declare(ticks=1.5);
declare(ticks=0);
declare(ticks=9223372036854775807);
declare(ticks=1e30);
declare(ticks="1e30");
declare(ticks="9999999999999999999999");
declare(ticks="12abc");
declare(ticks=0x7FFFFFFFFFFFFFFFF);
echo "compiled\n";
