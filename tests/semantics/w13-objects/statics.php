<?php
// REQ-018; T-035/036. Static cells and instance properties have distinct owners.
class W13StaticCells {public static int $p=1;public static int $u;public static ?int $n;protected static int $hidden=2;public int $instance=3;}
$tests['static-instance-read']=function(){$o=new W13StaticCells;return [$o->p,W13StaticCells::$p];};
$tests['static-instance-write']=function(){$o=new W13StaticCells;$o->p='dynamic';return [$o->p,W13StaticCells::$p];};
$tests['static-instance-reference']=function(){$o=new W13StaticCells;$r=&$o->p;$r='alias';return [$o->p,W13StaticCells::$p];};
$tests['static-instance-unset']=function(){$o=new W13StaticCells;unset($o->p);return W13StaticCells::$p;};
$tests['static-instance-isset']=function(){$o=new W13StaticCells;return [isset($o->p),isset(W13StaticCells::$p)];};
$tests['static-instance-protected']=function(){$o=new W13StaticCells;return attempt(fn()=>$o->hidden);};
$tests['static-reference-type']=function(){$r=&W13StaticCells::$p;$e=attempt(function()use(&$r){$r='bad';});return [$e,$r,W13StaticCells::$p];};
$tests['static-reference-uninitialized']=function(){return attempt(function(){$r=&W13StaticCells::$u;});};
$tests['static-reference-nullable']=function(){$r=&W13StaticCells::$n;return [$r,W13StaticCells::$n];};
$tests['static-unset']=function(){return [attempt(function(){unset(W13StaticCells::$p);}),attempt(function(){unset(W13StaticCells::$hidden);}),attempt(function(){unset(W13StaticCells::$missing);}),attempt(function(){unset(W13StaticCells::$instance);})];};
