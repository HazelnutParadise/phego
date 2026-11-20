<?php
function say($x) {
    echo "say ", $x, "\n";
    return $x;
}
echo "start\n";
no_such_function(...say([1]), a: say("never evaluated"));
