<?php
// finally_break_in_switch: a break inside a switch within try stays inside the switch; a return from a switch inside try runs finally
function pick($v)
{
    try {
        switch ($v) {
            case 1:
                echo "case 1\n";
                break;
            case 2:
                echo "case 2 then fallthrough\n";
            case 3:
                echo "case 3\n";
                break;
            default:
                echo "default\n";
        }
        echo "after switch for ", $v, "\n";
    } finally {
        echo "pick finally for ", $v, "\n";
    }
    return "picked " . $v;
}
$r = pick(1);
echo $r, "\n";
$r = pick(2);
echo $r, "\n";
$r = pick(9);
echo $r, "\n";

function returnFromSwitch($v)
{
    try {
        switch ($v) {
            case 'stop':
                return "returned from switch";
            default:
                echo "not stop\n";
        }
        return "after switch";
    } finally {
        echo "returnFromSwitch finally\n";
    }
}
$r = returnFromSwitch('stop');
echo $r, "\n";
$r = returnFromSwitch('go');
echo $r, "\n";

for ($i = 0; $i < 2; $i++) {
    try {
        switch ($i) {
            case 0:
                break;
        }
        echo "loop body ", $i, "\n";
    } finally {
        echo "loop finally ", $i, "\n";
    }
}
