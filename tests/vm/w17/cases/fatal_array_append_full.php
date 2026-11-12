<?php
$a = [9223372036854775807 => "max"];
echo "full\n";
$a[] = "overflow";
echo "never\n";
