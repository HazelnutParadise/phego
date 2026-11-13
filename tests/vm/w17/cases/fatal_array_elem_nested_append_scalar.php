<?php
$a = ["k" => 5];
echo "before\n";
$a["k"][] = 5;
echo "not reached\n";
