<?php
$s = "abc";
echo "before\n";
$s[0] .= "d";
echo "not reached\n";
