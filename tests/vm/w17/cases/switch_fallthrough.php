<?php
function sw($v)
{
    switch ($v) {
        case 1:
            echo "one ";
        case 2:
            echo "two ";
            break;
        default:
            echo "default ";
        case 3:
            echo "three ";
            break;
        case "abc":
            echo "abc ";
            break;
    }
    echo "|\n";
}
sw(1);
sw(2);
sw(3);
sw(4);
sw("1");
sw("1.0");
sw(true);
sw("abc");
sw(0);
sw(null);
switch ("") {
    case null:
        echo "empty matches null\n";
        break;
    case "":
        echo "never\n";
}
switch (true) {
    case 5 > 9:
        echo "never\n";
        break;
    case 5 > 2:
        echo "first true case\n";
        break;
}
switch (1) {
}
echo "end\n";
