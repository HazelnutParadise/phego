<?php
$i = 5;
echo "start\n";
$r = &$i[0];
echo "unreached\n";
