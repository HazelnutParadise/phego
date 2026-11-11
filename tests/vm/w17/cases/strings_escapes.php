<?php
echo 'single \'quoted\' \\ \n stays', "\n";
echo "tab[\t] newline-escape done\n";
echo "dollar \$x and backslash \\ and quote \"\n";
echo "hex \x41\x62 octal \101\060 unicode \u{48}\u{49}\n";
echo "unknown \q escape kept\n";
echo "vertical\v|escape\e|form\f|end\n";
echo 'multi
line single', "\n";
echo "\u{1F600}", "\n";
echo "nul[\0]byte\n";
