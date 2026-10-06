package deps

import (
	"strings"
	"testing"
)

func TestReadMap(t *testing.T) {
	src := `iwmap 4
"000_Global" flags expanded  active
"000_Global/No Comp" flags hidden ignore 
// entity 0
{
guid "{7C68F7A9}"
"classname" "worldspawn"
"skyboxmodel" "skybox_default_day"
// brush 0
{
 guid "{F8FC4ED4}"
 ( 0 0 -672 ) ( 5408 0 -672 ) ( 5408 32 -672 ) sky 16 16 2080 -2080 0 0 lightmap_gray 16384 16384 2080 -2080 0 0
 ( 4 32 1344 ) ( 5412 32 1344 ) ( 5412 0 1344 ) mtl_wall 16 16 2096 -2080 0 0 lightmap_gray 16384 16384 2096 -2080 0 0
}
// brush 2: in an ignored layer
 {
  layer "000_Global/No Comp"
  ( 0 0 0 ) ( 1 0 0 ) ( 1 1 0 ) mtl_hidden 16 16 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0
 }
// brush 1
 {
  guid "{9A94AB2F}"
  curve
  {
  toolFlags;
   mtl_rope
   lightmap_gray
   9 11 16 1
   (
	v 1175.2 -1555.5 129.2 t 0 0 1 1
   )
  }
 }
}
// entity 1
{
"classname" "misc_prefab"
"model" "_prefabs/zm/house.map"
}
// entity 2
{
layer "000_Global/No Comp/spawners"
"classname" "actor_zm_basic"
}
`
	ents, err := readMap(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 2 {
		t.Fatalf("%d entities (the one in an ignored layer is left out)", len(ents))
	}
	if got := strings.Join(ents[0].materials, ","); got != "sky,mtl_wall,mtl_rope" {
		t.Errorf("world materials: %s", got)
	}
	if ents[0].keys["skyboxmodel"] != "skybox_default_day" || ents[1].keys["model"] != "_prefabs/zm/house.map" {
		t.Errorf("keys: %v %v", ents[0].keys, ents[1].keys)
	}
}
