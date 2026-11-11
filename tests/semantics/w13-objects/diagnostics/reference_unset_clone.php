<?php
class H5 {public string $a='old';}
class HS5 {function __toString():string{unset($GLOBALS['r']);$GLOBALS['c']=clone$GLOBALS['o'];return 'new';}}
$GLOBALS['o']=new H5;$GLOBALS['r']=&$GLOBALS['o']->a;$GLOBALS['r']=new HS5;echo json_encode([$GLOBALS['o']->a,$GLOBALS['c']->a,array_key_exists('r',$GLOBALS)]),"\n";
