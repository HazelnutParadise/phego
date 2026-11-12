<?php
function inner($v)
{
    return 1.5 / $v;
}
function outer($v, $label)
{
    echo $label, "\n";
    return inner($v);
}
echo outer(0.0, "dividing"), "\n";
