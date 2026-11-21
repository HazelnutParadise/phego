<?php
namespace App\Models;

interface Model
{
    const TABLE = "none";
    public function table();
}

class User implements Model
{
    const TABLE = "users";
    public function table()
    {
        return static::TABLE;
    }
}

class Admin extends User
{
    const TABLE = "admins";
}

function describe(Model $m)
{
    return $m::class . " -> " . $m->table();
}

echo User::class, "\n";
echo \App\Models\User::class, "\n";
echo namespace\Admin::class, "\n";
echo Model::class, "\n";
echo describe(new User), "\n";
echo describe(new \App\Models\Admin), "\n";
$name = "App\\Models\\Admin";
$a = new $name;
echo "dynamic new: ", $a::class, " ", $a->table(), "\n";
$short = "Admin";
echo "unqualified string resolves globally: ", $short, "\n";
echo "instanceof qualified: ", ($a instanceof \App\Models\User ? "yes" : "no"), "\n";
echo "instanceof string: ", ($a instanceof $name ? "yes" : "no"), "\n";
echo "class name of unknown class constant: ", Missing\Thing::class, "\n";
echo "case kept as written: ", user::class, "\n";
echo __NAMESPACE__, "\n";
