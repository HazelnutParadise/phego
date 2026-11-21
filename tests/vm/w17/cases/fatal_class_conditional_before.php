<?php
echo "start\n";
$flag = true;
echo (new Maybe)->v, "\n";
if ($flag) {
    class Maybe
    {
        public $v = "declared";
    }
}
