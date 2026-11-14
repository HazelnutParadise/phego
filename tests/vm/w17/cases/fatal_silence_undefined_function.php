<?php
echo @$u1, "a\n";
echo @$u1 . $u2, "b\n";
function f()
{
    echo @$x;
    return 1;
}
echo f(), "c\n";
exit(@g());
