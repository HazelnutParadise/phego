<?php
function dup($v)
{
    return clone $v;
}
echo "cloning null\n";
dup(null);
