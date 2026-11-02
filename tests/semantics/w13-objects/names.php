<?php
// PHP folds ASCII letters, preserving non-ASCII identifier bytes.
class W13NameMethods {public function Ö(){return 'upper';}public function ö(){return 'lower';}}
class W13Ö {}
class W13ö {}
class W13NameTypes {public W13Ö $o;}
$tests['method-non-ascii']=function(){$o=new W13NameMethods;return [$o->Ö(),$o->ö()];};
$tests['class-non-ascii']=function(){return [get_class(new W13Ö),get_class(new W13ö)];};
$tests['property-non-ascii-type']=function(){$o=new W13NameTypes;return attempt(function()use($o){$o->o=new W13ö;});};
