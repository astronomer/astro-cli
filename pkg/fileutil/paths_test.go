package fileutil

func (s *Suite) TestGetWorkingDir() {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			name:    "basic case",
			want:    "fileutil",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := GetWorkingDir()
			if (err != nil) != tt.wantErr {
				s.Errorf(err, "GetWorkingDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			s.Contains(got, tt.want)
		})
	}
}

func (s *Suite) TestGetHomeDir() {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			name:    "basic case",
			want:    "/",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := GetHomeDir()
			if (err != nil) != tt.wantErr {
				s.Errorf(err, "GetHomeDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			s.Contains(got, tt.want)
		})
	}
}

func (s *Suite) TestIsEmptyDir() {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "basic case",
			args: args{path: "."},
			want: false,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(IsEmptyDir(tt.args.path), tt.want)
		})
	}
}
