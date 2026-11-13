<?php
$a = ["s" => "str"];
echo "before\n";
unset($a["s"][0]);
echo "not reached\n";
