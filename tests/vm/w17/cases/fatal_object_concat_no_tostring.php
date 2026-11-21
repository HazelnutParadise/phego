<?php
class Plain
{
}
function label($o)
{
    return "label: " . $o;
}
echo "labelling\n";
echo label(new Plain), "\n";
