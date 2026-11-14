<?php
function f()
{
    return @g();
}
function g()
{
    echo $u;
    return "g";
}
echo f(), "\n";
echo $v, "\n";
function h()
{
    @($x = $w);
    echo $y;
    return 1;
}
echo h(), "\n";
function loud()
{
    echo $loud_u;
    return "loud";
}
function quiet_caller()
{
    $r = @loud();
    echo $after_u;
    return $r;
}
echo quiet_caller(), "\n";
function returns_inside()
{
    echo @$a;
    return @(1 . $b);
}
echo returns_inside(), $c, "\n";
