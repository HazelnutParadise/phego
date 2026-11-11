<?php
function caller($v)
{
    echo "in caller\n";
    return Missing_Thing($v);
}
caller([1, 2]);
