<?php
// exception_destructor_timing: destruction of a user exception object around catch, unset, overwrite and function exit
class NoisyError extends Exception {
    public function __destruct() {
        echo "noisy destroyed: ", $this->getMessage(), "\n";
    }
}

echo "step1\n";
$x = new NoisyError("a");
$x = null;
echo "step2\n";

try {
    throw new NoisyError("b");
} catch (NoisyError $c) {
    echo "caught b\n";
}
unset($c);
echo "step3\n";

try {
    throw new NoisyError("c");
} catch (NoisyError $d) {
    echo "caught c\n";
}
try {
    throw new NoisyError("d");
} catch (NoisyError $d) {
    echo "caught d over c\n";
}
echo "step4\n";

function local_catch() {
    try {
        throw new NoisyError("e");
    } catch (NoisyError $l) {
        echo "in local_catch\n";
    }
    echo "local_catch end\n";
}
local_catch();
echo "after local_catch\n";

$keep = new NoisyError("f");
echo "end of script\n";
