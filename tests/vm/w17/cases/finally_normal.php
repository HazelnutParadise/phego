<?php
// finally_normal: try/finally and try/catch/finally with no exception: order of the blocks
function plain()
{
    echo "plain: try\n";
    try {
        echo "plain: body\n";
    } finally {
        echo "plain: finally\n";
    }
    echo "plain: after\n";
}
plain();

function withCatch()
{
    try {
        echo "catch: try\n";
    } catch (Exception $e) {
        echo "catch: never\n";
    } finally {
        echo "catch: finally\n";
    }
    echo "catch: after\n";
}
withCatch();

function valueThrough()
{
    try {
        $x = "value";
    } finally {
        echo "value finally runs\n";
    }
    return $x;
}
$r = valueThrough();
echo "returned: ", $r, "\n";

for ($i = 1; $i <= 2; $i++) {
    try {
        echo "loop try ", $i, "\n";
    } finally {
        echo "loop finally ", $i, "\n";
    }
}

$v = 1;
try {
    $v = 2;
} finally {
    $v = $v + 10;
}
echo "v after finally: ", $v, "\n";
echo "top-level done\n";
