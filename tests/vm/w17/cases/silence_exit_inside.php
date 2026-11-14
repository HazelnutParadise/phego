<?php
function f()
{
    echo $u;
    @g();
    echo $v;
}
function g()
{
    echo $w;
    exit("bye\n");
}
f();
