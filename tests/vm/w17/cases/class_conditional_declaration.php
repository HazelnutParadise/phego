<?php
$mode = "b";

if ($mode === "a") {
    class Impl
    {
        public function which()
        {
            return "A implementation";
        }
    }
} else {
    class Impl
    {
        public function which()
        {
            return "B implementation";
        }
    }
}
echo (new Impl)->which(), "\n";

function declare_inside()
{
    class FromFunction
    {
        public $v = "declared when the function ran";
    }
    echo "declared\n";
}
declare_inside();
echo (new FromFunction)->v, "\n";

$flag = true;
if ($flag) {
    class Only
    {
        const X = "only";
    }
}
echo Only::X, "\n";

$n = 0;
while ($n < 1) {
    class InLoop
    {
        const N = "loop class";
    }
    $n++;
}
echo InLoop::N, "\n";
